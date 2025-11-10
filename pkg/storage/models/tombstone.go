package models

import (
	"fmt"
	"time"
)

// Tombstone represents a deleted object marker
type Tombstone struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // OBJECT#{object_id}
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // TOMBSTONE

	// GSI keys for querying tombstones by actor and type
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"GSI1PK"` // ACTOR#{actor_id}#TOMBSTONES
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"GSI1SK"` // DELETED#{timestamp}

	// GSI for querying tombstones by type
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsI2PK" json:"GSI2PK"` // TOMBSTONE#{former_type}
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsI2SK" json:"GSI2SK"` // DELETED#{timestamp}

	// Core fields from legacy
	ID         string    `dynamorm:"attr:id" json:"id"`                          // Original object ID
	Type       string    `dynamorm:"attr:type" json:"type"`                      // Always "Tombstone"
	FormerType string    `dynamorm:"attr:formerType" json:"formerType"`          // Original object type
	Deleted    time.Time `dynamorm:"attr:deleted" json:"deleted"`                // When it was deleted
	DeletedBy  string    `dynamorm:"attr:deletedBy" json:"deletedBy"`            // Actor who deleted it
	Summary    string    `dynamorm:"attr:summary" json:"summary,omitempty"`      // Optional deletion reason
	CreatedAt  time.Time `dynamorm:"attr:createdAt" json:"CreatedAt"`            // When the tombstone was created

	// TTL field for automatic cleanup after 30 days
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl"`
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
	if err := t.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return nil
}

// UpdateKeys updates GSI keys for the tombstone
func (t *Tombstone) UpdateKeys() error {
	// Validate required fields
	if t.ID == "" {
		return fmt.Errorf("ID is required")
	}

	// Set primary keys
	t.PK = fmt.Sprintf(KeyPatternObject, t.ID)
	t.SK = "TOMBSTONE"

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
	return nil
}

// GetPK returns the primary key for BaseRepository interface
func (t *Tombstone) GetPK() string {
	return t.PK
}

// GetSK returns the sort key for BaseRepository interface
func (t *Tombstone) GetSK() string {
	return t.SK
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
