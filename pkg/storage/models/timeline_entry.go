package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// TimelineEntry represents an entry in a user's timeline stored in DynamoDB
type TimelineEntry struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // TIMELINE#{type}#{id}
	SK string `theorydb:"sk,attr:SK" json:"-"` // {timestamp}#{entryID}

	// GSI for public timeline queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"` // TIMELINE#PUBLIC#{local/federated}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"` // {timestamp}#{entryID}

	// Entry data
	TimelineType string    `theorydb:"attr:timelineType" json:"timeline_type"` // HOME, PUBLIC, LIST, DIRECT
	TimelineID   string    `theorydb:"attr:timelineID" json:"timeline_id"`     // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST
	EntryID      string    `theorydb:"attr:entryID" json:"entry_id"`           // Unique ID for this entry (usually timestamp + post ID)
	PostID       string    `theorydb:"attr:postID" json:"post_id"`             // The actual post/object ID
	ActorID      string    `theorydb:"attr:actorID" json:"actor_id"`           // Who created the post
	ActorHandle  string    `theorydb:"attr:actorHandle" json:"actor_handle"`   // Actor's handle for quick display
	Content      string    `theorydb:"attr:content" json:"content"`            // First 500 chars for preview
	ContentType  string    `theorydb:"attr:contentType" json:"content_type"`   // Note, Article, etc.
	HasMedia     bool      `theorydb:"attr:hasMedia" json:"has_media"`         // Quick flag for media
	IsReply      bool      `theorydb:"attr:isReply" json:"is_reply"`           // Is this a reply?
	InReplyTo    string    `theorydb:"attr:inReplyTo" json:"in_reply_to"`      // ID of post being replied to
	IsBoost      bool      `theorydb:"attr:isBoost" json:"is_boost"`           // Is this a boost/announce?
	BoostedBy    string    `theorydb:"attr:boostedBy" json:"boosted_by"`       // Who boosted it (if applicable)
	Visibility   string    `theorydb:"attr:visibility" json:"visibility"`      // public, unlisted, private, direct
	Language     string    `theorydb:"attr:language" json:"language"`          // Language code
	Sensitive    bool      `theorydb:"attr:sensitive" json:"sensitive"`        // Content warning flag
	SpoilerText  string    `theorydb:"attr:spoilerText" json:"spoiler_text"`   // Content warning text
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"created_at"`       // When the post was created
	TimelineAt   time.Time `theorydb:"attr:timelineAt" json:"timeline_at"`     // When it was added to timeline (for sorting)
	ExpiresAt    time.Time `theorydb:"attr:expiresAt" json:"expires_at"`       // TTL for auto-deletion

	// TTL for DynamoDB auto-deletion
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	if err := common.ValidateRequiredParam("e.EntryID", e.EntryID); err != nil {
		e.EntryID = fmt.Sprintf("%d#%s", e.TimelineAt.UnixNano(), e.PostID)
	}
}

// IsExpired checks if the timeline entry has expired
func (e *TimelineEntry) IsExpired() bool {
	return !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)
}

// TableName returns the DynamoDB table backing TimelineEntry.
func (TimelineEntry) TableName() string {
	return MainTableName
}
