package models

import (
	"fmt"
	"time"
)

// ReviewerStats represents statistics about a moderation reviewer
type ReviewerStats struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // REVIEWER#{reviewerID}
	SK string `theorydb:"sk,attr:SK" json:"-"` // STATS

	// Attributes from interface
	ReviewerID        string         `theorydb:"attr:reviewerID" json:"reviewer_id"`
	TotalReviews      int            `theorydb:"attr:totalReviews" json:"total_reviews"`
	AccurateReviews   int            `theorydb:"attr:accurateReviews" json:"accurate_reviews"`
	AccuracyRate      float64        `theorydb:"attr:accuracyRate" json:"accuracy_rate"`
	LastReviewAt      time.Time      `theorydb:"attr:lastReviewAt" json:"last_review_at"`
	TrustScore        float64        `theorydb:"attr:trustScore" json:"trust_score"`
	JoinedAt          time.Time      `theorydb:"attr:joinedAt" json:"joined_at"`
	ReviewsByCategory map[string]int `theorydb:"attr:reviewsByCategory" json:"reviews_by_category"`

	// Additional statistics
	ConsecutiveAccurate   int                `theorydb:"attr:consecutiveAccurate" json:"consecutive_accurate"`   // Current streak
	MaxStreak             int                `theorydb:"attr:maxStreak" json:"max_streak"`                       // Best streak
	RecentAccuracy        float64            `theorydb:"attr:recentAccuracy" json:"recent_accuracy"`             // Last 100 reviews
	ResponseTimeAvg       float64            `theorydb:"attr:responseTimeAvg" json:"response_time_avg"`          // Average response time in seconds
	DisagreementRate      float64            `theorydb:"attr:disagreementRate" json:"disagreement_rate"`         // Rate of disagreeing with consensus
	SpecializationScores  map[string]float64 `theorydb:"attr:specializationScores" json:"specialization_scores"` // Accuracy by category
	LastTrainingCompleted *time.Time         `theorydb:"attr:lastTrainingCompleted" json:"last_training_completed,omitempty"`
	BadgesEarned          []string           `theorydb:"attr:badgesEarned" json:"badges_earned,omitempty"`
	UpdatedAt             time.Time          `theorydb:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys updates the partition and sort keys
func (r *ReviewerStats) UpdateKeys() {
	r.PK = fmt.Sprintf("REVIEWER#%s", r.ReviewerID)
	r.SK = SKStats
}

// NewReviewerStats creates new reviewer statistics
func NewReviewerStats(reviewerID string) *ReviewerStats {
	stats := &ReviewerStats{
		ReviewerID:           reviewerID,
		JoinedAt:             time.Now().UTC(),
		LastReviewAt:         time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
		ReviewsByCategory:    make(map[string]int),
		SpecializationScores: make(map[string]float64),
		BadgesEarned:         []string{},
		TrustScore:           50.0, // Start at neutral trust
	}
	stats.UpdateKeys()
	return stats
}

// GetReviewerStatsKey returns the key for retrieving reviewer stats
func GetReviewerStatsKey(reviewerID string) (pk, sk string) {
	return fmt.Sprintf("REVIEWER#%s", reviewerID), "STATS"
}

// RecordReview updates stats after a review
func (r *ReviewerStats) RecordReview(category string, wasAccurate bool, responseTime float64) {
	r.TotalReviews++
	r.LastReviewAt = time.Now().UTC()
	r.UpdatedAt = time.Now().UTC()

	// Update category counts
	if r.ReviewsByCategory == nil {
		r.ReviewsByCategory = make(map[string]int)
	}
	r.ReviewsByCategory[category]++

	// Update accuracy
	if wasAccurate {
		r.AccurateReviews++
		r.ConsecutiveAccurate++
		if r.ConsecutiveAccurate > r.MaxStreak {
			r.MaxStreak = r.ConsecutiveAccurate
		}
	} else {
		r.ConsecutiveAccurate = 0
	}

	// Update accuracy rate
	r.CalculateAccuracyRate()

	// Update response time average
	if r.TotalReviews == 1 {
		r.ResponseTimeAvg = responseTime
	} else {
		r.ResponseTimeAvg = ((r.ResponseTimeAvg * float64(r.TotalReviews-1)) + responseTime) / float64(r.TotalReviews)
	}

	// Update trust score
	r.CalculateTrustScore()
}

// CalculateAccuracyRate calculates the overall accuracy rate
func (r *ReviewerStats) CalculateAccuracyRate() {
	if r.TotalReviews > 0 {
		r.AccuracyRate = (float64(r.AccurateReviews) / float64(r.TotalReviews)) * 100
	} else {
		r.AccuracyRate = 0
	}
}

// CalculateTrustScore calculates trust score based on multiple factors
func (r *ReviewerStats) CalculateTrustScore() {
	// Base score from accuracy (0-50 points)
	accuracyScore := r.AccuracyRate * 0.5

	// Experience bonus (0-20 points)
	experienceScore := 0.0
	if r.TotalReviews >= 1000 {
		experienceScore = 20.0
	} else if r.TotalReviews >= 500 {
		experienceScore = 15.0
	} else if r.TotalReviews >= 100 {
		experienceScore = 10.0
	} else if r.TotalReviews >= 50 {
		experienceScore = 5.0
	}

	// Consistency bonus (0-15 points)
	consistencyScore := 0.0
	if r.ConsecutiveAccurate >= 50 {
		consistencyScore = 15.0
	} else if r.ConsecutiveAccurate >= 25 {
		consistencyScore = 10.0
	} else if r.ConsecutiveAccurate >= 10 {
		consistencyScore = 5.0
	}

	// Speed bonus (0-10 points) - faster reviews get more points
	speedScore := 0.0
	if r.ResponseTimeAvg > 0 && r.ResponseTimeAvg < 30 {
		speedScore = 10.0
	} else if r.ResponseTimeAvg < 60 {
		speedScore = 7.0
	} else if r.ResponseTimeAvg < 120 {
		speedScore = 4.0
	}

	// Activity bonus (0-5 points) - recent activity
	activityScore := 0.0
	daysSinceLastReview := time.Since(r.LastReviewAt).Hours() / 24
	if daysSinceLastReview < 1 {
		activityScore = 5.0
	} else if daysSinceLastReview < 7 {
		activityScore = 3.0
	} else if daysSinceLastReview < 30 {
		activityScore = 1.0
	}

	r.TrustScore = accuracyScore + experienceScore + consistencyScore + speedScore + activityScore

	// Cap at 100
	if r.TrustScore > 100 {
		r.TrustScore = 100
	}
}

// UpdateSpecializationScore updates accuracy for a specific category
func (r *ReviewerStats) UpdateSpecializationScore(category string, recentAccuracy float64) {
	if r.SpecializationScores == nil {
		r.SpecializationScores = make(map[string]float64)
	}
	r.SpecializationScores[category] = recentAccuracy
}

// GetSpecialization returns the reviewer's best category
func (r *ReviewerStats) GetSpecialization() (category string, score float64) {
	for cat, acc := range r.SpecializationScores {
		if acc > score && r.ReviewsByCategory[cat] >= 50 { // Minimum 50 reviews
			category = cat
			score = acc
		}
	}
	return
}

// IsExperienced checks if reviewer has enough experience
func (r *ReviewerStats) IsExperienced() bool {
	return r.TotalReviews >= 100 && r.AccuracyRate >= 80
}

// IsTrusted checks if reviewer is highly trusted
func (r *ReviewerStats) IsTrusted() bool {
	return r.TrustScore >= 80
}

// NeedsTraining checks if reviewer needs additional training
func (r *ReviewerStats) NeedsTraining() bool {
	// Needs training if accuracy drops below 70% or hasn't trained in 90 days
	needsAccuracyTraining := r.AccuracyRate < 70 && r.TotalReviews >= 20

	needsRefresher := false
	if r.LastTrainingCompleted != nil {
		daysSinceTraining := time.Since(*r.LastTrainingCompleted).Hours() / 24
		needsRefresher = daysSinceTraining > 90
	}

	return needsAccuracyTraining || needsRefresher
}

// TableName returns the DynamoDB table backing ReviewerStats.
func (ReviewerStats) TableName() string {
	return MainTableName
}

// EarnBadge adds a badge if not already earned
func (r *ReviewerStats) EarnBadge(badge string) bool {
	for _, b := range r.BadgesEarned {
		if b == badge {
			return false // Already has badge
		}
	}
	r.BadgesEarned = append(r.BadgesEarned, badge)
	r.UpdatedAt = time.Now().UTC()
	return true
}
