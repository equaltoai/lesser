package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
)

// AIAnalysis represents an AI analysis result stored in DynamoDB
type AIAnalysis struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "AI#{object_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "ANALYSIS#{analysis_id}"

	// GSI4 for temporal queries (reusing cost tracking GSI)
	GSI4PK string `theorydb:"index:gsi4,pk,attr:gsi4PK" json:"gsi4_pk"` // Format: "AI#ANALYSIS#{date}"
	GSI4SK string `theorydb:"index:gsi4,sk,attr:gsi4SK" json:"gsi4_sk"` // Format: "{timestamp}"

	// Core fields
	ID         string    `theorydb:"attr:id" json:"id"`
	ObjectID   string    `theorydb:"attr:objectID" json:"object_id"`
	ObjectType string    `theorydb:"attr:objectType" json:"object_type"`
	AnalyzedAt time.Time `theorydb:"attr:analyzedAt" json:"analyzed_at"`
	Version    string    `theorydb:"attr:version" json:"version"`

	// Analysis results
	TextAnalysis  *ai.TextAnalysis  `theorydb:"attr:textAnalysis" json:"text_analysis,omitempty"`
	ImageAnalysis *ai.ImageAnalysis `theorydb:"attr:imageAnalysis" json:"image_analysis,omitempty"`
	AIDetection   *ai.AIDetection   `theorydb:"attr:aiDetection" json:"ai_detection,omitempty"`
	SpamAnalysis  *ai.SpamAnalysis  `theorydb:"attr:spamAnalysis" json:"spam_analysis,omitempty"`

	// Composite scores
	OverallRisk      float64 `theorydb:"attr:overallRisk" json:"overall_risk"`
	ModerationAction string  `theorydb:"attr:moderationAction" json:"moderation_action"`
	Confidence       float64 `theorydb:"attr:confidence" json:"confidence"`

	// DynamoDB metadata
	Type      string    `theorydb:"attr:type" json:"type"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Using object keys for updating
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "OBJECT#{object_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "OBJECT#{object_id}"

	// Queue metadata
	ForceAnalysis bool      `theorydb:"attr:forceAnalysis" json:"force_analysis"`
	UpdatedAt     time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
