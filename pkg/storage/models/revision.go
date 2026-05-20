package models

import (
	"fmt"
	"time"
)

// Revision represents a historical version of a published object
type Revision struct {
	// Primary keys: OBJECT#{object_id}#REVISION / VERSION#{version}
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// Core fields
	ID       string `theorydb:"attr:id" json:"id"` // Unique revision ID
	ObjectID string `theorydb:"attr:objectID" json:"object_id"`
	Version  int    `theorydb:"attr:version" json:"version"`

	// Snapshot of content at this version
	Content      string `theorydb:"attr:content" json:"content"`
	ContentHash  string `theorydb:"attr:contentHash" json:"content_hash"` // SHA256 for deduplication
	MetadataJSON string `theorydb:"attr:metadataJSON" json:"metadata_json"`

	// Change tracking
	ChangeSummary string `theorydb:"attr:changeSummary" json:"change_summary,omitempty"` // Optional commit message
	ChangedBy     string `theorydb:"attr:changedBy" json:"changed_by"`
	ChangeType    string `theorydb:"attr:changeType" json:"change_type"` // create, update, restore
	GeneratedBy   string `theorydb:"attr:generatedBy,omitempty" json:"generated_by,omitempty"`
	ReviewedBy    string `theorydb:"attr:reviewedBy,omitempty" json:"reviewed_by,omitempty"`
	PublishedBy   string `theorydb:"attr:publishedBy,omitempty" json:"published_by,omitempty"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
