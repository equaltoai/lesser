package models

import (
	"fmt"
	"time"
)

// Revision represents a historical version of a published object
type Revision struct {
	// Primary keys: OBJECT#{object_id}#REVISION / VERSION#{version}
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// Core fields
	ID       string `dynamorm:"attr:id" json:"id"` // Unique revision ID
	ObjectID string `dynamorm:"attr:objectID" json:"object_id"`
	Version  int    `dynamorm:"attr:version" json:"version"`

	// Snapshot of content at this version
	Content      string `dynamorm:"attr:content" json:"content"`
	ContentHash  string `dynamorm:"attr:contentHash" json:"content_hash"` // SHA256 for deduplication
	MetadataJSON string `dynamorm:"attr:metadataJSON" json:"metadata_json"`

	// Change tracking
	ChangeSummary string `dynamorm:"attr:changeSummary" json:"change_summary,omitempty"` // Optional commit message
	ChangedBy     string `dynamorm:"attr:changedBy" json:"changed_by"`
	ChangeType    string `dynamorm:"attr:changeType" json:"change_type"` // create, update, restore

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing Revision.
func (Revision) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Revision model
func (r *Revision) UpdateKeys() error {
	if r.ObjectID == "" {
		return fmt.Errorf("ObjectID is required")
	}

	r.PK = fmt.Sprintf("OBJECT#%s#REVISION", r.ObjectID)
	r.SK = fmt.Sprintf("VERSION#%08d", r.Version) // Zero-padded for sort order

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	r.UpdatedAt = r.CreatedAt

	return nil
}

// GetPK returns the partition key
func (r *Revision) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *Revision) GetSK() string {
	return r.SK
}
