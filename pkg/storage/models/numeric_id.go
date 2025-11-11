package models

import (
	"time"
)

// NumericIDMapping represents a mapping from numeric ID to username for Mastodon API compatibility
type NumericIDMapping struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "NUMERIC_ID#{numeric_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "METADATA"

	// Mapping data
	NumericID string    `dynamorm:"attr:numericID" json:"numeric_id"`
	Username  string    `dynamorm:"attr:username" json:"username"`
	ActorID   string    `dynamorm:"attr:actorID" json:"actor_id"`
	Type      string    `dynamorm:"attr:type" json:"type"` // "NumericIDMapping"
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table name for the NumericIDMapping model
func (NumericIDMapping) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (n *NumericIDMapping) BeforeCreate() error {
	n.PK = "NUMERIC_ID#" + n.NumericID
	n.SK = SKMetadata
	n.Type = "NumericIDMapping"
	n.CreatedAt = time.Now()
	return nil
}
