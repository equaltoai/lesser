package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
)

// AIAnalysis represents an AI analysis result stored in DynamoDB
type AIAnalysis struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "AI#{object_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "ANALYSIS#{analysis_id}"

	// GSI4 for temporal queries (reusing cost tracking GSI)
	GSI4PK string `dynamorm:"index:cost-date-index,pk,attr:gsI4PK" json:"gsi4_pk"` // Format: "AI#ANALYSIS#{date}"
	GSI4SK string `dynamorm:"index:cost-date-index,sk,attr:gsI4SK" json:"gsi4_sk"` // Format: "{timestamp}"

	// Core fields
	ID         string    `dynamorm:"attr:id" json:"id"`
	ObjectID   string    `dynamorm:"attr:objectID" json:"object_id"`
	ObjectType string    `dynamorm:"attr:objectType" json:"object_type"`
	AnalyzedAt time.Time `dynamorm:"attr:analyzedAt" json:"analyzed_at"`
	Version    string    `dynamorm:"attr:version" json:"version"`

	// Analysis results
	TextAnalysis  *ai.TextAnalysis  `dynamorm:"attr:textAnalysis" json:"text_analysis,omitempty"`
	ImageAnalysis *ai.ImageAnalysis `dynamorm:"attr:imageAnalysis" json:"image_analysis,omitempty"`
	AIDetection   *ai.AIDetection   `dynamorm:"attr:aiDetection" json:"ai_detection,omitempty"`
	SpamAnalysis  *ai.SpamAnalysis  `dynamorm:"attr:spamAnalysis" json:"spam_analysis,omitempty"`

	// Composite scores
	OverallRisk      float64 `dynamorm:"attr:overallRisk" json:"overall_risk"`
	ModerationAction string  `dynamorm:"attr:moderationAction" json:"moderation_action"`
	Confidence       float64 `dynamorm:"attr:confidence" json:"confidence"`

	// DynamoDB metadata
	Type      string    `dynamorm:"attr:type" json:"type"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys updates the GSI keys for the AI analysis
func (a *AIAnalysis) UpdateKeys() error {
	a.PK = "AI#" + a.ObjectID
	a.SK = "ANALYSIS#" + a.ID
	a.GSI4PK = "AI#ANALYSIS#" + a.AnalyzedAt.Format(common.DateFormat)
	a.GSI4SK = a.AnalyzedAt.Format(time.RFC3339Nano)
	a.Type = "AIAnalysis"
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (a *AIAnalysis) GetPK() string {
	return a.PK
}

// GetSK returns the sort key for BaseModel interface
func (a *AIAnalysis) GetSK() string {
	return a.SK
}

// BeforeCreate hook for DynamORM
func (a *AIAnalysis) BeforeCreate() error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	a.UpdatedAt = time.Now()
	return a.UpdateKeys()
}

// BeforeUpdate hook for DynamORM
func (a *AIAnalysis) BeforeUpdate() error {
	a.UpdatedAt = time.Now()
	return a.UpdateKeys()
}

// TableName returns the DynamoDB table backing AIAnalysis.
func (AIAnalysis) TableName() string {
	return MainTableName
}

// AIAnalysisQueue represents a queued object for AI analysis
type AIAnalysisQueue struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Using object keys for updating
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "OBJECT#{object_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "OBJECT#{object_id}"

	// Queue metadata
	ForceAnalysis bool      `dynamorm:"attr:forceAnalysis" json:"force_analysis"`
	UpdatedAt     time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys updates the keys for the queue entry
func (q *AIAnalysisQueue) UpdateKeys() error {
	// Keys are set externally since they depend on the object ID
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (q *AIAnalysisQueue) GetPK() string {
	return q.PK
}

// GetSK returns the sort key for BaseModel interface
func (q *AIAnalysisQueue) GetSK() string {
	return q.SK
}

// BeforeUpdate hook for DynamORM
func (q *AIAnalysisQueue) BeforeUpdate() error {
	q.UpdatedAt = time.Now()
	return q.UpdateKeys()
}

// TableName returns the DynamoDB table backing AIAnalysisQueue.
func (AIAnalysisQueue) TableName() string {
	return MainTableName
}
