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

	// GSI keys for querying tombstones by actor and type
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"` // ACTOR#{actor_id}#TOMBSTONES  
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK"` // DELETED#{timestamp}

	// GSI for querying tombstones by type
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"GSI2PK"` // TOMBSTONE#{former_type}
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"GSI2SK"` // DELETED#{timestamp}

	// Core fields from legacy
	ID         string    `json:"id"`                // Original object ID
	Type       string    `json:"type"`              // Always "Tombstone"
	FormerType string    `json:"formerType"`        // Original object type
	Deleted    time.Time `json:"deleted"`           // When it was deleted
	DeletedBy  string    `json:"deletedBy"`         // Actor who deleted it
	Summary    string    `json:"summary,omitempty"` // Optional deletion reason
	CreatedAt  time.Time `json:"CreatedAt"`         // When the tombstone was created
	
	// TTL field for automatic cleanup after 30 days
	TTL int64 `dynamorm:"ttl" json:"ttl"`
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

	// Set TTL to 30 days from now (for automatic cleanup)
	if t.TTL == 0 {
		t.TTL = time.Now().AddDate(0, 0, 30).Unix()
	}

	// Set keys
	t.PK = fmt.Sprintf(KeyPatternObject, t.ID)
	t.SK = "TOMBSTONE"

	// Update GSI keys
	t.UpdateKeys()

	return nil
}

// UpdateKeys updates GSI keys for the tombstone
func (t *Tombstone) UpdateKeys() {
	// GSI1: Query tombstones by actor
	if t.DeletedBy != "" {
		t.GSI1PK = fmt.Sprintf("ACTOR#%s#TOMBSTONES", t.DeletedBy)
		t.GSI1SK = fmt.Sprintf("DELETED#%d", t.Deleted.Unix())
	}

	// GSI2: Query tombstones by former type
	if t.FormerType != "" {
		t.GSI2PK = fmt.Sprintf("TOMBSTONE#%s", t.FormerType)
		t.GSI2SK = fmt.Sprintf("DELETED#%d", t.Deleted.Unix())
	}
}

// IsTombstone always returns true for tombstone objects
func (t *Tombstone) IsTombstone() bool {
	return true
}

// GetOriginalID returns the ID of the original object
func (t *Tombstone) GetOriginalID() string {
	return t.ID
}

// GetDeletedBy returns the actor who deleted the object
func (t *Tombstone) GetDeletedBy() string {
	return t.DeletedBy
}

// GetFormerType returns the type of the original object
func (t *Tombstone) GetFormerType() string {
	return t.FormerType
}

// ShouldCleanup returns true if the tombstone is past its TTL
func (t *Tombstone) ShouldCleanup() bool {
	return time.Now().Unix() > t.TTL
}
