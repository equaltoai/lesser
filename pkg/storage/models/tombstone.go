package models

import (
	"fmt"
	"time"
)

// Tombstone represents a deleted object marker
type Tombstone struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // OBJECT#{object_id}
	SK string `dynamorm:"sk" json:"SK"` // TOMBSTONE

	// Core fields from legacy
	ID         string    `json:"id"`                // Original object ID
	Type       string    `json:"type"`              // Always "Tombstone"
	FormerType string    `json:"formerType"`        // Original object type
	Deleted    time.Time `json:"deleted"`           // When it was deleted
	DeletedBy  string    `json:"deletedBy"`         // Actor who deleted it
	Summary    string    `json:"summary,omitempty"` // Optional deletion reason
	CreatedAt  time.Time `json:"CreatedAt"`         // When the tombstone was created
}

// TableName returns the DynamoDB table name
func (Tombstone) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the Tombstone for creation
func (t *Tombstone) BeforeCreate() error {
	// Set type
	t.Type = "Tombstone"

	// Set timestamps if not already set
	if t.Deleted.IsZero() {
		t.Deleted = time.Now()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	// Set keys
	t.PK = fmt.Sprintf(KeyPatternObject, t.ID)
	t.SK = "TOMBSTONE"

	return nil
}
