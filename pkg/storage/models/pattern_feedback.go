package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// PatternFeedback represents feedback on pattern matching results
type PatternFeedback struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // PATTERN#{patternID}
	SK string `theorydb:"sk,attr:SK" json:"-"` // FEEDBACK#{timestamp}#{feedbackID}

	// Attributes from interface
	WasMatch         bool `theorydb:"attr:wasMatch" json:"was_match"`
	WasFalsePositive bool `theorydb:"attr:wasFalsePositive" json:"was_false_positive"`

	// Additional attributes
	PatternID   string    `theorydb:"attr:patternID" json:"pattern_id"`
	FeedbackID  string    `theorydb:"attr:feedbackID" json:"feedback_id"`
	SubmittedBy string    `theorydb:"attr:submittedBy" json:"submitted_by"` // User or system that submitted feedback
	SubmittedAt time.Time `theorydb:"attr:submittedAt" json:"submitted_at"`
	ContentID   string    `theorydb:"attr:contentID" json:"content_id"`     // ID of content that was evaluated
	ContentType string    `theorydb:"attr:contentType" json:"content_type"` // Type of content (status, user, etc)
	PatternType string    `theorydb:"attr:patternType" json:"pattern_type"` // spam, abuse, etc
	Confidence  float64   `theorydb:"attr:confidence" json:"confidence"`    // Original confidence score
	Notes       string    `theorydb:"attr:notes" json:"notes,omitempty"`    // Additional feedback notes
	TTL         int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`    // 90 days retention
}

// UpdateKeys updates the partition and sort keys
func (p *PatternFeedback) UpdateKeys() {
	p.PK = fmt.Sprintf("PATTERN#%s", p.PatternID)
	p.SK = fmt.Sprintf("FEEDBACK#%s#%s", p.SubmittedAt.Format(time.RFC3339Nano), p.FeedbackID)

	// Set TTL to 90 days from submission
	p.TTL = p.SubmittedAt.AddDate(0, 3, 0).Unix()
}

// NewPatternFeedback creates a new pattern feedback entry
func NewPatternFeedback(patternID, contentID, submittedBy string) *PatternFeedback {
	feedback := &PatternFeedback{
		PatternID:   patternID,
		FeedbackID:  uuid.New().String(),
		ContentID:   contentID,
		SubmittedBy: submittedBy,
		SubmittedAt: time.Now().UTC(),
	}
	feedback.UpdateKeys()
	return feedback
}

// GetPatternFeedbackKey returns the key for retrieving specific feedback
func GetPatternFeedbackKey(patternID, timestamp, feedbackID string) (pk, sk string) {
	return fmt.Sprintf("PATTERN#%s", patternID),
		fmt.Sprintf("FEEDBACK#%s#%s", timestamp, feedbackID)
}

// GetPatternFeedbackKeys returns keys for querying all feedback for a pattern
func GetPatternFeedbackKeys(patternID string) (pk, skPrefix string) {
	return fmt.Sprintf("PATTERN#%s", patternID), "FEEDBACK#"
}

// GetPatternFeedbackRangeKeys returns keys for querying feedback in a time range
func GetPatternFeedbackRangeKeys(patternID string, startTime, endTime time.Time) (pk, skStart, skEnd string) {
	pk = fmt.Sprintf("PATTERN#%s", patternID)
	skStart = fmt.Sprintf("FEEDBACK#%s", startTime.Format(time.RFC3339Nano))
	skEnd = fmt.Sprintf("FEEDBACK#%s", endTime.Format(time.RFC3339Nano))
	return
}

// IsCorrect returns true if the pattern matched correctly
func (p *PatternFeedback) IsCorrect() bool {
	// Pattern was correct if:
	// 1. It matched and wasn't a false positive, OR
	// 2. It didn't match and this was the right decision
	return (p.WasMatch && !p.WasFalsePositive) || (!p.WasMatch && p.WasFalsePositive)
}

// GetFeedbackType returns a string describing the type of feedback
func (p *PatternFeedback) GetFeedbackType() string {
	switch {
	case p.WasMatch && !p.WasFalsePositive:
		return "true_positive"
	case p.WasMatch && p.WasFalsePositive:
		return "false_positive"
	case !p.WasMatch && p.WasFalsePositive:
		return "false_negative"
	default:
		return "true_negative"
	}
}

// CalculatePatternAccuracy calculates accuracy from a slice of feedback
func CalculatePatternAccuracy(feedbacks []*PatternFeedback) float64 {
	if err := common.ValidateSliceNotEmpty("feedbacks", feedbacks); err != nil {
		return 0
	}

	correct := 0
	for _, f := range feedbacks {
		if f.IsCorrect() {
			correct++
		}
	}

	return float64(correct) / float64(len(feedbacks)) * 100
}

// CalculatePatternMetrics calculates detailed metrics from feedback
func CalculatePatternMetrics(feedbacks []*PatternFeedback) map[string]interface{} {
	metrics := map[string]interface{}{
		"total_feedback":  len(feedbacks),
		"true_positives":  0,
		"false_positives": 0,
		"true_negatives":  0,
		"false_negatives": 0,
		"accuracy":        0.0,
		"precision":       0.0,
		"recall":          0.0,
	}

	if err := common.ValidateSliceNotEmpty("feedbacks", feedbacks); err != nil {
		return metrics
	}

	tp, fp, tn, fn := 0, 0, 0, 0

	for _, f := range feedbacks {
		switch f.GetFeedbackType() {
		case "true_positive":
			tp++
		case "false_positive":
			fp++
		case "true_negative":
			tn++
		case "false_negative":
			fn++
		}
	}

	metrics["true_positives"] = tp
	metrics["false_positives"] = fp
	metrics["true_negatives"] = tn
	metrics["false_negatives"] = fn

	// Calculate accuracy
	if total := tp + fp + tn + fn; total > 0 {
		metrics["accuracy"] = float64(tp+tn) / float64(total) * 100
	}

	// Calculate precision (how many selected items are relevant)
	if tp+fp > 0 {
		metrics["precision"] = float64(tp) / float64(tp+fp) * 100
	}

	// Calculate recall (how many relevant items are selected)
	if tp+fn > 0 {
		metrics["recall"] = float64(tp) / float64(tp+fn) * 100
	}

	return metrics
}

// TableName returns the DynamoDB table backing PatternFeedback.
func (PatternFeedback) TableName() string {
	return MainTableName
}
