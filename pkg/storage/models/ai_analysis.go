package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
)

// AIAnalysis represents an AI analysis result stored in DynamoDB
type AIAnalysis struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "AI#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "ANALYSIS#{analysis_id}"

	// GSI4 for temporal queries (reusing cost tracking GSI)
	GSI4PK string `dynamorm:"index:cost-date-index,pk" json:"gsi4_pk"` // Format: "AI#ANALYSIS#{date}"
	GSI4SK string `dynamorm:"index:cost-date-index,sk" json:"gsi4_sk"` // Format: "{timestamp}"

	// Core fields
	ID         string    `json:"id"`
	ObjectID   string    `json:"object_id"`
	ObjectType string    `json:"object_type"`
	AnalyzedAt time.Time `json:"analyzed_at"`
	Version    string    `json:"version"`

	// Analysis results
	TextAnalysis  *ai.TextAnalysis  `json:"text_analysis,omitempty"`
	ImageAnalysis *ai.ImageAnalysis `json:"image_analysis,omitempty"`
	AIDetection   *ai.AIDetection   `json:"ai_detection,omitempty"`
	SpamAnalysis  *ai.SpamAnalysis  `json:"spam_analysis,omitempty"`

	// Composite scores
	OverallRisk      float64 `json:"overall_risk"`
	ModerationAction string  `json:"moderation_action"`
	Confidence       float64 `json:"confidence"`

	// DynamoDB metadata
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateKeys updates the GSI keys for the AI analysis
func (a *AIAnalysis) UpdateKeys() {
	a.PK = "AI#" + a.ObjectID
	a.SK = "ANALYSIS#" + a.ID
	a.GSI4PK = "AI#ANALYSIS#" + a.AnalyzedAt.Format(common.DateFormat)
	a.GSI4SK = a.AnalyzedAt.Format(time.RFC3339Nano)
	a.Type = "AIAnalysis"
}

// BeforeCreate hook for DynamORM
func (a *AIAnalysis) BeforeCreate() error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	a.UpdatedAt = time.Now()
	a.UpdateKeys()
	return nil
}

// BeforeUpdate hook for DynamORM
func (a *AIAnalysis) BeforeUpdate() error {
	a.UpdatedAt = time.Now()
	a.UpdateKeys()
	return nil
}

// AIAnalysisQueue represents a queued object for AI analysis
type AIAnalysisQueue struct {
	// Using object keys for updating
	PK string `dynamorm:"pk" json:"pk"` // Format: "OBJECT#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "OBJECT#{object_id}"

	// Queue metadata
	ForceAnalysis bool      `json:"force_analysis"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// UpdateKeys updates the keys for the queue entry
func (q *AIAnalysisQueue) UpdateKeys() {
	// Keys are set externally since they depend on the object ID
}

// BeforeUpdate hook for DynamORM
func (q *AIAnalysisQueue) BeforeUpdate() error {
	q.UpdatedAt = time.Now()
	return nil
}
