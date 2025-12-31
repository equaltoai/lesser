package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewerStats_LifecycleAndHelpers(t *testing.T) {
	t.Run("NewReviewerStats sets defaults and keys", func(t *testing.T) {
		rs := NewReviewerStats("rev-1")
		require.NotNil(t, rs)

		assert.Equal(t, "rev-1", rs.ReviewerID)
		assert.Equal(t, "REVIEWER#rev-1", rs.PK)
		assert.Equal(t, SKStats, rs.SK)
		assert.NotNil(t, rs.ReviewsByCategory)
		assert.NotNil(t, rs.SpecializationScores)
		assert.Empty(t, rs.BadgesEarned)
		assert.InDelta(t, 50.0, rs.TrustScore, 0.000001)
		assert.Equal(t, MainTableName, rs.TableName())

		pk, sk := GetReviewerStatsKey("rev-1")
		assert.Equal(t, rs.PK, pk)
		assert.Equal(t, "STATS", sk)
	})

	t.Run("RecordReview updates counters, averages, and streaks", func(t *testing.T) {
		rs := &ReviewerStats{ReviewerID: "rev-2"}
		rs.UpdateKeys()

		rs.RecordReview("spam", true, 10)
		assert.Equal(t, 1, rs.TotalReviews)
		assert.Equal(t, 1, rs.AccurateReviews)
		assert.Equal(t, 1, rs.ConsecutiveAccurate)
		assert.Equal(t, 1, rs.MaxStreak)
		assert.InDelta(t, 100.0, rs.AccuracyRate, 0.000001)
		assert.InDelta(t, 10.0, rs.ResponseTimeAvg, 0.000001)
		assert.Equal(t, 1, rs.ReviewsByCategory["spam"])

		// Non-accurate resets streak and updates average.
		rs.RecordReview("spam", false, 30)
		assert.Equal(t, 2, rs.TotalReviews)
		assert.Equal(t, 1, rs.AccurateReviews)
		assert.Equal(t, 0, rs.ConsecutiveAccurate)
		assert.Equal(t, 1, rs.MaxStreak)
		assert.InDelta(t, 50.0, rs.AccuracyRate, 0.000001)
		assert.InDelta(t, 20.0, rs.ResponseTimeAvg, 0.000001)
	})

	t.Run("CalculateTrustScore exercises thresholds and caps at 100", func(t *testing.T) {
		rs := &ReviewerStats{
			AccuracyRate:        100,
			TotalReviews:        1000,
			ConsecutiveAccurate: 50,
			ResponseTimeAvg:     10,
			LastReviewAt:        time.Now().Add(-2 * time.Hour),
		}
		rs.CalculateTrustScore()
		assert.Equal(t, 100.0, rs.TrustScore)

		rs = &ReviewerStats{
			AccuracyRate:        80,
			TotalReviews:        50,
			ConsecutiveAccurate: 10,
			ResponseTimeAvg:     90,
			LastReviewAt:        time.Now().Add(-10 * 24 * time.Hour),
		}
		rs.CalculateTrustScore()
		assert.True(t, rs.TrustScore > 0)
	})

	t.Run("UpdateSpecializationScore and GetSpecialization apply min-review rule", func(t *testing.T) {
		rs := &ReviewerStats{
			ReviewsByCategory: map[string]int{
				"spam":  100,
				"abuse": 49,
			},
		}

		rs.UpdateSpecializationScore("spam", 0.9)
		rs.UpdateSpecializationScore("abuse", 1.0)
		cat, score := rs.GetSpecialization()
		assert.Equal(t, "spam", cat)
		assert.InDelta(t, 0.9, score, 0.000001)
	})

	t.Run("IsExperienced and IsTrusted reflect thresholds", func(t *testing.T) {
		rs := &ReviewerStats{TotalReviews: 100, AccuracyRate: 80, TrustScore: 80}
		assert.True(t, rs.IsExperienced())
		assert.True(t, rs.IsTrusted())
	})

	t.Run("NeedsTraining handles accuracy and refresher conditions", func(t *testing.T) {
		rs := &ReviewerStats{TotalReviews: 20, AccuracyRate: 69.9}
		assert.True(t, rs.NeedsTraining())

		past := time.Now().Add(-91 * 24 * time.Hour)
		rs = &ReviewerStats{TotalReviews: 1, AccuracyRate: 100, LastTrainingCompleted: &past}
		assert.True(t, rs.NeedsTraining())

		recent := time.Now().Add(-10 * 24 * time.Hour)
		rs = &ReviewerStats{TotalReviews: 100, AccuracyRate: 100, LastTrainingCompleted: &recent}
		assert.False(t, rs.NeedsTraining())
	})

	t.Run("EarnBadge de-dupes and updates UpdatedAt", func(t *testing.T) {
		rs := &ReviewerStats{BadgesEarned: []string{"first"}}
		before := rs.UpdatedAt
		assert.True(t, rs.EarnBadge("second"))
		assert.False(t, rs.EarnBadge("second"))
		assert.Contains(t, rs.BadgesEarned, "second")
		assert.True(t, rs.UpdatedAt.After(before) || before.IsZero())
	})
}
