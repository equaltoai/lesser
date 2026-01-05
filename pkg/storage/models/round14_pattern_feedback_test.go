package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternFeedback_KeysTypesAndAggregations(t *testing.T) {
	t.Run("NewPatternFeedback and UpdateKeys set keys and TTL", func(t *testing.T) {
		pf := NewPatternFeedback("p1", "c1", "system")
		require.NotNil(t, pf)
		assert.Equal(t, "PATTERN#p1", pf.PK)
		assert.Contains(t, pf.SK, "FEEDBACK#")
		assert.NotEmpty(t, pf.FeedbackID)
		assert.True(t, pf.TTL > 0)
		assert.Equal(t, MainTableName, (PatternFeedback{}).TableName())
	})

	t.Run("Key helpers", func(t *testing.T) {
		pk, sk := GetPatternFeedbackKey("p1", "ts", "id")
		assert.Equal(t, "PATTERN#p1", pk)
		assert.Equal(t, "FEEDBACK#ts#id", sk)

		pk, prefix := GetPatternFeedbackKeys("p1")
		assert.Equal(t, "PATTERN#p1", pk)
		assert.Equal(t, "FEEDBACK#", prefix)

		start := time.Unix(1700000000, 0).UTC()
		end := start.Add(time.Hour)
		pk, skStart, skEnd := GetPatternFeedbackRangeKeys("p1", start, end)
		assert.Equal(t, "PATTERN#p1", pk)
		assert.Contains(t, skStart, "FEEDBACK#")
		assert.Contains(t, skEnd, "FEEDBACK#")
	})

	t.Run("IsCorrect and GetFeedbackType cover truth table", func(t *testing.T) {
		pf := &PatternFeedback{WasMatch: true, WasFalsePositive: false}
		assert.True(t, pf.IsCorrect())
		assert.Equal(t, "true_positive", pf.GetFeedbackType())

		pf = &PatternFeedback{WasMatch: true, WasFalsePositive: true}
		assert.False(t, pf.IsCorrect())
		assert.Equal(t, "false_positive", pf.GetFeedbackType())

		pf = &PatternFeedback{WasMatch: false, WasFalsePositive: true}
		assert.True(t, pf.IsCorrect())
		assert.Equal(t, "false_negative", pf.GetFeedbackType())

		pf = &PatternFeedback{WasMatch: false, WasFalsePositive: false}
		assert.False(t, pf.IsCorrect())
		assert.Equal(t, "true_negative", pf.GetFeedbackType())
	})

	t.Run("CalculatePatternAccuracy returns percent", func(t *testing.T) {
		assert.Equal(t, 0.0, CalculatePatternAccuracy(nil))

		feedbacks := []*PatternFeedback{
			{WasMatch: true, WasFalsePositive: false},  // correct
			{WasMatch: true, WasFalsePositive: true},   // incorrect
			{WasMatch: false, WasFalsePositive: true},  // correct
			{WasMatch: false, WasFalsePositive: false}, // incorrect
		}
		assert.InDelta(t, 50.0, CalculatePatternAccuracy(feedbacks), 0.000001)
	})

	t.Run("CalculatePatternMetrics computes counts and rates", func(t *testing.T) {
		empty := CalculatePatternMetrics(nil)
		assert.Equal(t, 0, empty["total_feedback"])

		feedbacks := []*PatternFeedback{
			{WasMatch: true, WasFalsePositive: false},  // TP
			{WasMatch: true, WasFalsePositive: true},   // FP
			{WasMatch: false, WasFalsePositive: true},  // FN (as modeled)
			{WasMatch: false, WasFalsePositive: false}, // TN
		}

		metrics := CalculatePatternMetrics(feedbacks)
		assert.Equal(t, 4, metrics["total_feedback"])
		assert.Equal(t, 1, metrics["true_positives"])
		assert.Equal(t, 1, metrics["false_positives"])
		assert.Equal(t, 1, metrics["true_negatives"])
		assert.Equal(t, 1, metrics["false_negatives"])

		assert.InDelta(t, 50.0, metrics["accuracy"].(float64), 0.000001)
		assert.InDelta(t, 50.0, metrics["precision"].(float64), 0.000001)
		assert.InDelta(t, 50.0, metrics["recall"].(float64), 0.000001)
	})
}

