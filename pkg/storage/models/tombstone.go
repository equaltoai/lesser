package models

import (
	"fmt"
	"time"
)

// Tombstone represents a deleted object marker
type Tombstone struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"PK"` // OBJECT#{object_id}
	SK string `theorydb:"sk,attr:SK" json:"SK"` // TOMBSTONE

	// GSI keys for querying tombstones by actor and type
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK"` // ACTOR#{actor_id}#TOMBSTONES
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK"` // DELETED#{timestamp}

	// GSI for querying tombstones by type
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2PK"` // TOMBSTONE#{former_type}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2SK"` // DELETED#{timestamp}

	// Core fields from legacy
	ID           string    `theorydb:"attr:id" json:"id"`                             // Original object ID
	Type         string    `theorydb:"attr:type" json:"type"`                         // Always "Tombstone"
	FormerType   string    `theorydb:"attr:formerType" json:"formerType"`             // Original object type
	Deleted      time.Time `theorydb:"attr:deleted" json:"deleted"`                   // When it was deleted
	DeletedBy    string    `theorydb:"attr:deletedBy" json:"deletedBy"`               // Actor who deleted it
	AttributedTo string    `theorydb:"attr:attributedTo,omitempty" json:"attributedTo,omitempty"` // Original object's author actor ID
	Summary      string    `theorydb:"attr:summary" json:"summary,omitempty"`         // Optional deletion reason
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`               // When the tombstone was created

	// TTL field for automatic cleanup after 30 days
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
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
