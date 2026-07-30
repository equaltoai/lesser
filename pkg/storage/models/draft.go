package models

import (
	"fmt"
	"strings"
	"time"
)

// Draft represents an unpublished content draft
type Draft struct {
	// Primary keys: USER#{author_id}#DRAFT / ID#{draft_id}
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI1: Object drafts - OBJECT#{object_id}#DRAFT / TIME#{updated_at}
	// Allows finding all drafts for a specific published object
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	// GSI4: Scheduled publishing + status index - DRAFT#STATUS#{status} / TIME#{timestamp}#AUTHOR#{author_id}#ID#{draft_id}
	// Allows finding drafts by status, and enables scheduled publishing workers to query due drafts efficiently.
	GSI4PK string `theorydb:"index:gsi4,pk,attr:gsi4PK,omitempty"`
	GSI4SK string `theorydb:"index:gsi4,sk,attr:gsi4SK,omitempty"`

	// Core fields
	ID       string  `theorydb:"attr:id" json:"id"`
	AuthorID string  `theorydb:"attr:authorID" json:"author_id"`
	ObjectID *string `theorydb:"attr:objectID" json:"object_id,omitempty"` // nil = new, set = editing existing

	// Content
	ContentType   string `theorydb:"attr:contentType" json:"content_type"` // Note, Article
	Title         string `theorydb:"attr:title" json:"title,omitempty"`    // For Article
	Slug          string `theorydb:"attr:slug" json:"slug,omitempty"`
	Content       string `theorydb:"attr:content" json:"content"`
	ContentFormat string `theorydb:"attr:contentFormat" json:"content_format"` // html, markdown

	// Draft state
	Status      string     `theorydb:"attr:status" json:"status"` // draft, scheduled, publishing, failed
	ScheduledAt *time.Time `theorydb:"attr:scheduledAt" json:"scheduled_at,omitempty"`

	// Metadata snapshot (full object metadata for preview)
	MetadataJSON string `theorydb:"attr:metadataJSON" json:"metadata_json,omitempty"`

	// Authoring attribution
	GeneratedBy  string `theorydb:"attr:generatedBy,omitempty" json:"generated_by,omitempty"`
	ReviewedBy   string `theorydb:"attr:reviewedBy,omitempty" json:"reviewed_by,omitempty"`
	ReviewStatus string `theorydb:"attr:reviewStatus,omitempty" json:"review_status,omitempty"`
	EditorNotes  string `theorydb:"attr:editorNotes,omitempty" json:"editor_notes,omitempty"`

	// Autosave tracking
	AutosaveVersion int       `theorydb:"attr:autosaveVersion" json:"autosave_version"`
	LastSavedAt     time.Time `theorydb:"attr:lastSavedAt" json:"last_saved_at"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
