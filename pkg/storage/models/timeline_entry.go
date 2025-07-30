package models

import (
	"fmt"
	"time"
)

// TimelineEntry represents an entry in a user's timeline stored in DynamoDB
type TimelineEntry struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // TIMELINE#{type}#{id}
	SK string `dynamorm:"sk" json:"-"` // {timestamp}#{entryID}

	// GSI for public timeline queries
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // TIMELINE#PUBLIC#{local/federated}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // {timestamp}#{entryID}

	// Entry data
	TimelineType string    `json:"timeline_type"` // HOME, PUBLIC, LIST, DIRECT
	TimelineID   string    `json:"timeline_id"`   // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST
	EntryID      string    `json:"entry_id"`      // Unique ID for this entry (usually timestamp + post ID)
	PostID       string    `json:"post_id"`       // The actual post/object ID
	ActorID      string    `json:"actor_id"`      // Who created the post
	ActorHandle  string    `json:"actor_handle"`  // Actor's handle for quick display
	Content      string    `json:"content"`       // First 500 chars for preview
	ContentType  string    `json:"content_type"`  // Note, Article, etc.
	HasMedia     bool      `json:"has_media"`     // Quick flag for media
	IsReply      bool      `json:"is_reply"`      // Is this a reply?
	InReplyTo    string    `json:"in_reply_to"`   // ID of post being replied to
	IsBoost      bool      `json:"is_boost"`      // Is this a boost/announce?
	BoostedBy    string    `json:"boosted_by"`    // Who boosted it (if applicable)
	Visibility   string    `json:"visibility"`    // public, unlisted, private, direct
	Language     string    `json:"language"`      // Language code
	Sensitive    bool      `json:"sensitive"`     // Content warning flag
	SpoilerText  string    `json:"spoiler_text"`  // Content warning text
	CreatedAt    time.Time `json:"created_at"`    // When the post was created
	TimelineAt   time.Time `json:"timeline_at"`   // When it was added to timeline (for sorting)
	ExpiresAt    time.Time `json:"expires_at"`    // TTL for auto-deletion

	// TTL for DynamoDB auto-deletion
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the timeline entry data
func (e *TimelineEntry) UpdateKeys() {
	// Primary key
	e.PK = fmt.Sprintf("TIMELINE#%s#%s", e.TimelineType, e.TimelineID)
	e.SK = fmt.Sprintf("%d#%s", e.TimelineAt.Unix(), e.EntryID)

	// GSI for public timeline
	if e.TimelineType == "PUBLIC" {
		e.GSI1PK = fmt.Sprintf("TIMELINE#PUBLIC#%s", e.TimelineID) // LOCAL or FEDERATED
		e.GSI1SK = e.SK
	} else {
		e.GSI1PK = ""
		e.GSI1SK = ""
	}

	// Set TTL if ExpiresAt is set
	if !e.ExpiresAt.IsZero() {
		e.TTL = e.ExpiresAt.Unix()
	}
}

// SetEntryID generates a unique entry ID if not already set
func (e *TimelineEntry) SetEntryID() {
	if e.EntryID == "" {
		e.EntryID = fmt.Sprintf("%d#%s", e.TimelineAt.UnixNano(), e.PostID)
	}
}

// IsExpired checks if the timeline entry has expired
func (e *TimelineEntry) IsExpired() bool {
	return !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)
}