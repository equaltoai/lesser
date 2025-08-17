// Package reputation provides actor reputation calculation algorithms based on activity history and trust metrics.
package reputation

import (
	"context"
	"math"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// Calculator computes reputation scores for actors
type Calculator struct {
	store       core.RepositoryStorage
	logger      *zap.Logger
	instanceURL string
}

// NewCalculator creates a new reputation calculator
func NewCalculator(store core.RepositoryStorage, instanceURL string, logger *zap.Logger) *Calculator {
	return &Calculator{
		store:       store,
		logger:      logger,
		instanceURL: instanceURL,
	}
}

// Calculate computes a reputation score for an actor
func (c *Calculator) Calculate(_ context.Context, input *CalculationInput) (*Reputation, error) {
	rep := &Reputation{
		ActorID:      input.ActorID,
		InstanceURL:  c.instanceURL, // Use configured instance URL
		CalculatedAt: time.Now(),
		Version:      "1.0",

		// Evidence
		TotalPosts:     input.PostCount,
		TotalFollowers: input.FollowerCount,
		AccountAge:     int(time.Since(input.AccountCreated).Hours() / 24),
		VouchCount:     len(input.VouchesReceived),
	}

	// Calculate individual scores
	rep.TrustScore = c.calculateTrustScore(input)
	rep.ActivityScore = c.calculateActivityScore(input)
	rep.ModerationScore = c.calculateModerationScore(input)
	rep.CommunityScore = c.calculateCommunityScore(input)

	// Total score is sum of all scores (max 1000)
	rep.TotalScore = rep.TrustScore + rep.ActivityScore + rep.ModerationScore + rep.CommunityScore

	c.logger.Info("Calculated reputation",
		zap.String("actor", input.ActorID),
		zap.Int("total_score", rep.TotalScore),
		zap.Int("trust", rep.TrustScore),
		zap.Int("activity", rep.ActivityScore),
		zap.Int("moderation", rep.ModerationScore),
		zap.Int("community", rep.CommunityScore))

	return rep, nil
}

// calculateTrustScore computes trust score based on trust graph (0-250)
func (c *Calculator) calculateTrustScore(input *CalculationInput) int {
	if err := common.ValidateSliceNotEmpty("input.TrustRelationships", input.TrustRelationships); err != nil {
		return 0
	}

	var totalTrust float64
	var weightedTrust float64
	recentRelationships := 0

	for _, rel := range input.TrustRelationships {
		// Only count incoming trust
		if rel.ToActor != input.ActorID {
			continue
		}

		// Weight by recency
		age := time.Since(rel.UpdatedAt)
		recencyWeight := 1.0
		if age > 30*24*time.Hour {
			recencyWeight = 0.5
		} else if age > 90*24*time.Hour {
			recencyWeight = 0.25
		}

		totalTrust += rel.TrustScore
		weightedTrust += rel.TrustScore * recencyWeight

		if age < 30*24*time.Hour {
			recentRelationships++
		}
	}

	// Base score on weighted average
	avgTrust := 0.0
	if len(input.TrustRelationships) > 0 {
		avgTrust = weightedTrust / float64(len(input.TrustRelationships))
	}

	// Scale to 0-200 based on average trust
	baseScore := int(avgTrust * 200)

	// Bonus for number of recent trusting relationships (up to 50)
	diversityBonus := int(math.Min(float64(recentRelationships*10), 50))

	score := baseScore + diversityBonus
	if score > 250 {
		score = 250
	}

	return score
}

// calculateActivityScore computes activity score (0-250)
func (c *Calculator) calculateActivityScore(input *CalculationInput) int {
	score := 0

	// Account age (up to 50 points)
	ageDays := int(time.Since(input.AccountCreated).Hours() / 24)
	ageScore := int(math.Min(float64(ageDays)/365*50, 50))
	score += ageScore

	// Post frequency (up to 100 points)
	postsPerDay := 0.0
	if ageDays > 0 {
		postsPerDay = float64(input.PostCount) / float64(ageDays)
	}

	// Ideal is 1-5 posts per day
	postScore := 0
	if postsPerDay >= 0.1 && postsPerDay <= 10 {
		if postsPerDay <= 5 {
			postScore = int(postsPerDay * 20) // Max 100 at 5 posts/day
		} else {
			postScore = 100 - int((postsPerDay-5)*10) // Penalty for over-posting
		}
	}
	score += postScore

	// Follower count (up to 50 points)
	followerScore := int(math.Min(math.Log10(float64(input.FollowerCount+1))*25, 50))
	score += followerScore

	// Recency bonus (up to 50 points)
	daysSinceActive := int(time.Since(input.LastActive).Hours() / 24)
	recencyScore := 0
	if daysSinceActive < 7 {
		recencyScore = 50
	} else if daysSinceActive < 30 {
		recencyScore = 30
	} else if daysSinceActive < 90 {
		recencyScore = 10
	}
	score += recencyScore

	if score > 250 {
		score = 250
	}

	return score
}

// calculateModerationScore computes moderation score (0-250)
func (c *Calculator) calculateModerationScore(input *CalculationInput) int {
	// Start with full score
	score := 250

	// Analyze moderation history
	reportsUpheld := 0
	reportsDismissed := 0
	suspensions := 0

	for _, event := range input.ModerationHistory {
		switch event.Type {
		case "report":
			switch event.Outcome {
			case OutcomeUpheld:
				reportsUpheld++
				// Deduct based on severity
				score -= event.Severity * 10
			case OutcomeDismissed:
				reportsDismissed++
				// Bonus for false reports
				score += 5
			}
		case "suspension":
			suspensions++
			score -= 100
		}
	}

	// Account age factor - newer accounts lose more points
	ageDays := int(time.Since(input.AccountCreated).Hours() / 24)
	if ageDays < 30 && reportsUpheld > 0 {
		score -= 50 // Extra penalty for new accounts with violations
	}

	// Ensure score stays in bounds
	if score < 0 {
		score = 0
	} else if score > 250 {
		score = 250
	}

	return score
}

// calculateCommunityScore computes community contribution score (0-250)
func (c *Calculator) calculateCommunityScore(input *CalculationInput) int {
	score := 0

	// Vouches received (up to 100 points)
	vouchScore := 0
	for _, vouch := range input.VouchesReceived {
		if vouch.Active && !vouch.Revoked && time.Now().Before(vouch.ExpiresAt) {
			// Weight by voucher reputation and confidence
			contribution := float64(vouch.VoucherReputation) / 1000.0 * vouch.Confidence * 20
			vouchScore += int(contribution)
		}
	}
	if vouchScore > 100 {
		vouchScore = 100
	}
	score += vouchScore

	// Vouches given that weren't revoked (up to 50 points)
	goodVouches := 0
	for _, vouch := range input.VouchesGiven {
		if vouch.Active && !vouch.Revoked {
			goodVouches++
		}
	}
	vouchGivenScore := int(math.Min(float64(goodVouches*10), 50))
	score += vouchGivenScore

	// Community notes (up to 50 points)
	noteScore := int(math.Min(float64(input.CommunityNotes*5), 50))
	score += noteScore

	// Helpful votes (up to 50 points)
	helpfulScore := int(math.Min(float64(input.HelpfulVotes), 50))
	score += helpfulScore

	if score > 250 {
		score = 250
	}

	return score
}

// GetBoostFromVouches calculates initial reputation boost from vouches
func (c *Calculator) GetBoostFromVouches(vouches []Vouch) int {
	boost := 0

	for _, vouch := range vouches {
		if vouch.Active && !vouch.Revoked && time.Now().Before(vouch.ExpiresAt) {
			// Calculate boost: voucher reputation * confidence / 10
			contribution := float64(vouch.VoucherReputation) * vouch.Confidence / 10.0
			boost += int(contribution)
		}
	}

	// Cap at 200 points
	if boost > 200 {
		boost = 200
	}

	return boost
}
