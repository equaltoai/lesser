package models

import (
	"time"
)

// NumericIDMapping represents a mapping from numeric ID to username for Mastodon API compatibility
type NumericIDMapping struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "NUMERIC_ID#{numeric_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "METADATA"

	// Mapping data
	NumericID string    `json:"numeric_id"`
	Username  string    `json:"username"`
	ActorID   string    `json:"actor_id"`
	Type      string    `json:"type"` // "NumericIDMapping"
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name for the NumericIDMapping model
func (NumericIDMapping) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (n *NumericIDMapping) BeforeCreate() error {
	n.PK = "NUMERIC_ID#" + n.NumericID
	n.SK = "METADATA"
	n.Type = "NumericIDMapping"
	n.CreatedAt = time.Now()
	return nil
}