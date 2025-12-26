package models

import (
	"fmt"
	"strings"
	"time"
)

// Draft represents an unpublished content draft
type Draft struct {
	// Primary keys: USER#{author_id}#DRAFT / ID#{draft_id}
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// GSI1: Object drafts - OBJECT#{object_id}#DRAFT / TIME#{updated_at}
	// Allows finding all drafts for a specific published object
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`

	// GSI4: Scheduled publishing + status index - DRAFT#STATUS#{status} / TIME#{timestamp}#AUTHOR#{author_id}#ID#{draft_id}
	// Allows finding drafts by status, and enables scheduled publishing workers to query due drafts efficiently.
	GSI4PK string `dynamorm:"index:gsi4,pk,attr:gsi4PK"`
	GSI4SK string `dynamorm:"index:gsi4,sk,attr:gsi4SK"`

	// Core fields
	ID       string  `dynamorm:"attr:id" json:"id"`
	AuthorID string  `dynamorm:"attr:authorID" json:"author_id"`
	ObjectID *string `dynamorm:"attr:objectID" json:"object_id,omitempty"` // nil = new, set = editing existing

	// Content
	ContentType   string `dynamorm:"attr:contentType" json:"content_type"` // Note, Article
	Title         string `dynamorm:"attr:title" json:"title,omitempty"`    // For Article
	Slug          string `dynamorm:"attr:slug" json:"slug,omitempty"`
	Content       string `dynamorm:"attr:content" json:"content"`
	ContentFormat string `dynamorm:"attr:contentFormat" json:"content_format"` // html, markdown

	// Draft state
	Status      string     `dynamorm:"attr:status" json:"status"` // draft, scheduled, publishing, failed
	ScheduledAt *time.Time `dynamorm:"attr:scheduledAt" json:"scheduled_at,omitempty"`

	// Metadata snapshot (full object metadata for preview)
	MetadataJSON string `dynamorm:"attr:metadataJSON" json:"metadata_json,omitempty"`

	// Autosave tracking
	AutosaveVersion int       `dynamorm:"attr:autosaveVersion" json:"autosave_version"`
	LastSavedAt     time.Time `dynamorm:"attr:lastSavedAt" json:"last_saved_at"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing Draft.
func (Draft) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Draft model
func (d *Draft) UpdateKeys() error {
	if d.AuthorID == "" {
		return fmt.Errorf("AuthorID is required")
	}
	if d.ID == "" {
		return fmt.Errorf("ID is required")
	}

	d.PK = fmt.Sprintf("USER#%s#DRAFT", d.AuthorID)
	d.SK = fmt.Sprintf("ID#%s", d.ID)

	if d.ObjectID != nil && *d.ObjectID != "" {
		d.GSI1PK = fmt.Sprintf("OBJECT#%s#DRAFT", *d.ObjectID)
	} else {
		d.GSI1PK = fmt.Sprintf("USER#%s#NEWDRAFT", d.AuthorID)
	}

	// Ensure timestamps
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = time.Now()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}

	d.GSI1SK = fmt.Sprintf("TIME#%s", d.UpdatedAt.Format(time.RFC3339Nano))

	status := strings.ToLower(strings.TrimSpace(d.Status))
	if status == "" {
		status = "draft"
	}

	timestamp := d.UpdatedAt.UTC()
	if status == "scheduled" {
		if d.ScheduledAt != nil && !d.ScheduledAt.IsZero() {
			timestamp = d.ScheduledAt.UTC()
		} else {
			timestamp = time.Now().UTC()
		}
	}

	d.GSI4PK = fmt.Sprintf("DRAFT#STATUS#%s", status)
	d.GSI4SK = fmt.Sprintf("TIME#%s#AUTHOR#%s#ID#%s", timestamp.Format(time.RFC3339Nano), d.AuthorID, d.ID)

	return nil
}

// GetPK returns the partition key
func (d *Draft) GetPK() string {
	return d.PK
}

// GetSK returns the sort key
func (d *Draft) GetSK() string {
	return d.SK
}
