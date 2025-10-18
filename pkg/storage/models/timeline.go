package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Timeline represents an entry in a user's timeline stored in DynamoDB using DynamORM
type Timeline struct {
	// Primary key - using composite key for timeline entries
	PK string `dynamorm:"pk" json:"pk"` // Format: "timeline#{timeline_type}#{timeline_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "{timeline_at_timestamp}#{entry_id}"

	// GSI1 - Post timeline (all timeline entries for a specific post)
	GSI1PK string `dynamorm:"index:post-timeline-index,pk" json:"gsi1_pk"` // Format: "POST#{post_id}"
	GSI1SK string `dynamorm:"index:post-timeline-index,sk" json:"gsi1_sk"` // Format: "{timeline_at_timestamp}#{entry_id}"

	// GSI2 - Actor timeline (all timeline entries by an actor)
	GSI2PK string `dynamorm:"index:actor-timeline-index,pk" json:"gsi2_pk"` // Format: "ACTOR#{actor_id}"
	GSI2SK string `dynamorm:"index:actor-timeline-index,sk" json:"gsi2_sk"` // Format: "{timeline_at_timestamp}#{entry_id}"

	// GSI3 - Visibility timeline (entries by visibility level)
	GSI3PK string `dynamorm:"index:visibility-timeline-index,pk" json:"gsi3_pk,omitempty"` // Format: "VISIBILITY#{visibility}"
	GSI3SK string `dynamorm:"index:visibility-timeline-index,sk" json:"gsi3_sk,omitempty"` // Format: "{timeline_at_timestamp}#{entry_id}"

	// GSI4 - Language timeline (entries by language)
	GSI4PK string `dynamorm:"index:language-timeline-index,pk" json:"gsi4_pk,omitempty"` // Format: "LANGUAGE#{language}"
	GSI4SK string `dynamorm:"index:language-timeline-index,sk" json:"gsi4_sk,omitempty"` // Format: "{timeline_at_timestamp}#{entry_id}"

	// Core timeline data
	TimelineType string `json:"timeline_type"` // HOME, PUBLIC, LIST, DIRECT, HASHTAG
	TimelineID   string `json:"timeline_id"`   // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST, hashtag for HASHTAG
	EntryID      string `json:"entry_id"`      // Unique ID for this entry (usually timestamp + post ID)
	PostID       string `json:"post_id"`       // The actual post/object ID
	ActorID      string `json:"actor_id"`      // Who created the post
	ActorHandle  string `json:"actor_handle"`  // Actor's handle for quick display

	// Content preview for performance
	Content     string `json:"content"`      // First 500 chars for preview
	ContentType string `json:"content_type"` // Note, Article, etc.

	// Quick flags for filtering
	HasMedia    bool   `json:"has_media"`    // Quick flag for media
	IsReply     bool   `json:"is_reply"`     // Is this a reply?
	InReplyTo   string `json:"in_reply_to"`  // ID of post being replied to
	IsBoost     bool   `json:"is_boost"`     // Is this a boost/announce?
	BoostedBy   string `json:"boosted_by"`   // Who boosted it (if applicable)
	Visibility  string `json:"visibility"`   // public, unlisted, private, direct
	Language    string `json:"language"`     // Language code
	Sensitive   bool   `json:"sensitive"`    // Content warning flag
	SpoilerText string `json:"spoiler_text"` // Content warning text

	// Timestamps
	CreatedAt  time.Time `json:"created_at"`                   // When the post was created
	TimelineAt time.Time `json:"timeline_at"`                  // When it was added to timeline (for sorting)
	TTL        int64     `dynamorm:"ttl" json:"ttl,omitempty"` // DynamoDB TTL (Unix timestamp)

	// DynamORM metadata
	ModifiedAt time.Time `dynamorm:"updated_at" json:"modified_at"`
	Version    int       `dynamorm:"version" json:"version"`
}

// TableName returns the DynamoDB table name for the Timeline model
func (Timeline) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (t *Timeline) BeforeCreate() error {
	now := time.Now()
	t.ModifiedAt = now

	// Set timeline time if not already set
	if t.TimelineAt.IsZero() {
		t.TimelineAt = now
	}

	// Set created time if not already set
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}

	// Generate entry ID if not set
	if err := common.ValidateRequiredParam("t.EntryID", t.EntryID); err != nil {
		t.EntryID = fmt.Sprintf("%d_%s", t.TimelineAt.Unix(), t.PostID)
	}

	// Set up primary key - match legacy pattern
	t.PK = fmt.Sprintf("TIMELINE#%s#%s", t.TimelineType, t.TimelineID)
	// Use reverse timestamp for newest-first ordering like legacy
	reverseTimestamp := 9999999999 - t.TimelineAt.Unix()
	t.SK = fmt.Sprintf("%010d#%s", reverseTimestamp, t.PostID)

	// Set up GSI keys
	t.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (t *Timeline) BeforeUpdate() error {
	t.ModifiedAt = time.Now()

	// Update GSI keys in case indexed fields changed
	t.setupGSIKeys()

	return nil
}

// setupGSIKeys configures all GSI partition and sort keys
func (t *Timeline) setupGSIKeys() {
	entryID := t.EntryID
	timestamp := t.TimelineAt.Unix()
	reverseTimestamp := 9999999999 - timestamp
	timestampStr := fmt.Sprintf("%010d", reverseTimestamp)

	// GSI1 - Used for public timeline in legacy (TIMELINE#PUBLIC#LOCAL/FEDERATED)
	if t.TimelineType == TimelinePublic {
		t.GSI1PK = fmt.Sprintf("TIMELINE#PUBLIC#%s", t.TimelineID) // LOCAL or FEDERATED
		t.GSI1SK = fmt.Sprintf("%s#%s", timestampStr, t.PostID)
	} else if t.PostID != "" {
		// Also used for post timeline
		t.GSI1PK = "POST#" + t.PostID
		t.GSI1SK = fmt.Sprintf("%s#%s", timestampStr, entryID)
	}

	// GSI2 - Actor timeline
	if t.ActorID != "" {
		t.GSI2PK = "ACTOR#" + t.ActorID
		t.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, entryID)
	}

	// GSI3 - Visibility timeline (only for public/unlisted content)
	if t.Visibility == "public" || t.Visibility == "unlisted" {
		t.GSI3PK = "VISIBILITY#" + t.Visibility
		t.GSI3SK = fmt.Sprintf("%s#%s", timestampStr, entryID)
	} else {
		t.GSI3PK = ""
		t.GSI3SK = ""
	}

	// GSI4 - Language timeline (only if language is specified)
	if t.Language != "" {
		t.GSI4PK = "LANGUAGE#" + t.Language
		t.GSI4SK = fmt.Sprintf("%s#%s", timestampStr, entryID)
	} else {
		t.GSI4PK = ""
		t.GSI4SK = ""
	}

}

// IsExpired returns true if the timeline entry has expired
func (t *Timeline) IsExpired() bool {
	return t.TTL > 0 && time.Now().Unix() > t.TTL
}

// SetTTL sets the TTL for this timeline entry
func (t *Timeline) SetTTL(expiresAt time.Time) {
	if !expiresAt.IsZero() {
		t.TTL = expiresAt.Unix()
	}
}

// IsPublic returns true if the timeline entry is publicly visible
func (t *Timeline) IsPublic() bool {
	return t.Visibility == "public"
}

// IsUnlisted returns true if the timeline entry is unlisted
func (t *Timeline) IsUnlisted() bool {
	return t.Visibility == "unlisted"
}

// IsPrivate returns true if the timeline entry is private
func (t *Timeline) IsPrivate() bool {
	return t.Visibility == "private"
}

// IsDirect returns true if the timeline entry is a direct message
func (t *Timeline) IsDirect() bool {
	return t.Visibility == "direct"
}

// IsHomeTimeline returns true if this is a home timeline entry
func (t *Timeline) IsHomeTimeline() bool {
	return t.TimelineType == "HOME"
}

// IsPublicTimeline returns true if this is a public timeline entry
func (t *Timeline) IsPublicTimeline() bool {
	return t.TimelineType == TimelinePublic
}

// IsListTimeline returns true if this is a list timeline entry
func (t *Timeline) IsListTimeline() bool {
	return t.TimelineType == "LIST"
}

// IsDirectTimeline returns true if this is a direct timeline entry
func (t *Timeline) IsDirectTimeline() bool {
	return t.TimelineType == "DIRECT"
}

// IsHashtagTimeline returns true if this is a hashtag timeline entry
func (t *Timeline) IsHashtagTimeline() bool {
	return t.TimelineType == "HASHTAG"
}

// GetTimelineKey returns the timeline key for this entry
func (t *Timeline) GetTimelineKey() string {
	return fmt.Sprintf("%s#%s", t.TimelineType, t.TimelineID)
}

// GetSortKey returns the sort key for this entry
func (t *Timeline) GetSortKey() string {
	// Use reverse timestamp like legacy
	reverseTimestamp := 9999999999 - t.TimelineAt.Unix()
	return fmt.Sprintf("%010d#%s", reverseTimestamp, t.EntryID)
}

// UpdateKeys updates the GSI keys for this timeline entry (required by DynamORM)
func (t *Timeline) UpdateKeys() error {
	t.setupGSIKeys()
	return nil
}

// GetPK returns the partition key for this timeline entry (required by BaseModel interface)
func (t *Timeline) GetPK() string {
	return t.PK
}

// GetSK returns the sort key for this timeline entry (required by BaseModel interface)
func (t *Timeline) GetSK() string {
	return t.SK
}
